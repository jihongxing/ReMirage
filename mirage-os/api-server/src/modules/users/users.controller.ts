import {
  Controller,
  Get,
  Patch,
  Param,
  Query,
  Body,
  UseGuards,
} from '@nestjs/common';
import { UsersService } from './users.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { RolesGuard } from '../auth/roles.guard';
import { OwnerGuard } from '../auth/owner.guard';
import { Permissions } from '../auth/permissions.decorator';
import { Permission } from '../auth/rbac-matrix';
import { PaginationDto } from '../../common/pagination.dto';

@Controller('users')
@UseGuards(JwtAuthGuard, RolesGuard, OwnerGuard)
export class UsersController {
  constructor(private usersService: UsersService) {}

  @Get()
  @Permissions(Permission.USER_READ)
  findAll(@Query() pagination: PaginationDto) {
    return this.usersService.findAll(pagination.page, pagination.limit);
  }

  @Get(':id')
  @Permissions(Permission.USER_READ)
  findOne(@Param('id') id: string) {
    return this.usersService.findOne(id);
  }

  @Patch(':id/pubkey')
  @Permissions(Permission.USER_WRITE)
  bindPubkey(@Param('id') id: string, @Body('pubkey') pubkey: string) {
    return this.usersService.bindPubkey(id, pubkey);
  }

  @Patch(':id/deactivate')
  @Permissions(Permission.USER_WRITE)
  deactivate(@Param('id') id: string) {
    return this.usersService.deactivate(id);
  }
}
