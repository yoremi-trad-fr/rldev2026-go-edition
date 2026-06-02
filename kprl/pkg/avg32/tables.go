package avg32

var textWinNames = map[byte]string{
	0x01: "text_window_hide",
	0x02: "text_window_hide_effect",
	0x03: "text_window_hide_redraw",
	0x04: "text_window_mouse_wait",
	0x05: "text_window_clear",
}

var jumpSceneNames = map[byte]string{
	0x01: "jump_scene",
	0x02: "call_scene",
}

var formattedTextNames = map[byte]string{
	0x01: "draw_value",
	0x02: "draw_value_zero_padded",
	0x03: "draw_text_pointer",
	0x11: "draw_value_unknown1",
	0x13: "draw_value_unknown2",
}

var fadeNames = map[byte]string{
	0x01: "fade",
	0x02: "fade_timed",
	0x03: "fade_color",
	0x04: "fade_timed_color",
	0x10: "fill_screen",
	0x11: "fill_screen_color",
}

var waitNames = map[byte]string{
	0x01: "wait",
	0x02: "wait_mouse",
	0x03: "wait_set_base",
	0x04: "wait_from_base",
	0x05: "wait_from_base_mouse",
	0x06: "wait_set_base_value",
	0x10: "wait_0x10",
	0x11: "wait_0x11",
	0x12: "wait_0x12",
	0x13: "wait_0x13",
}

var graphicsNames = map[byte]string{
	0x01: "grp_load",
	0x02: "grp_load_effect",
	0x03: "grp_load2",
	0x04: "grp_load_effect2",
	0x05: "grp_load3",
	0x06: "grp_load_effect3",
	0x08: "grp_unknown1",
	0x09: "grp_load_to_buffer",
	0x10: "grp_load_to_buffer2",
	0x11: "grp_load_caching",
	0x13: "grp_cmd_0x13",
	0x22: "grp_load_composite",
	0x24: "grp_load_composite_indexed",
	0x30: "macro_buffer_clear",
	0x31: "macro_buffer_delete",
	0x32: "macro_buffer_read",
	0x33: "macro_buffer_set",
	0x50: "backup_screen_copy",
	0x52: "backup_screen_display",
	0x54: "grp_load_to_buffer3",
}

var soundNames = map[byte]string{
	0x01: "bgm_loop",
	0x02: "bgm_wait",
	0x03: "bgm_once",
	0x05: "bgm_fadein_loop",
	0x06: "bgm_fadein_wait",
	0x07: "bgm_fadein_once",
	0x10: "bgm_fadeout",
	0x11: "bgm_stop",
	0x12: "bgm_rewind",
	0x16: "bgm_unknown1",
	0x20: "koe_play_wait",
	0x21: "koe_play",
	0x22: "koe_play2",
	0x30: "wav_play",
	0x31: "wav_play2",
	0x32: "wav_loop",
	0x33: "wav_loop2",
	0x34: "wav_play_wait",
	0x35: "wav_play_wait2",
	0x36: "wav_stop",
	0x37: "wav_stop2",
	0x38: "wav_stop3",
	0x39: "wav_unknown_0x39",
	0x40: "se_play",
	0x50: "movie_play",
	0x51: "movie_loop",
	0x52: "movie_wait",
	0x53: "movie_wait_cancelable",
	0x54: "movie_wait2",
	0x55: "movie_wait_cancelable2",
	0x60: "sound_unknown1",
}

var choiceNames = map[byte]string{
	0x01: "choice",
	0x02: "choice2",
	0x04: "load_menu",
}

var stringNames = map[byte]string{
	0x01: "strcpy_literal",
	0x02: "strlen",
	0x03: "strcmp",
	0x04: "strcat",
	0x05: "strcpy",
	0x06: "itoa",
	0x07: "han_to_zen",
	0x08: "atoi",
}

var setMultiNames = map[byte]string{
	0x01: "set_multi_value",
	0x02: "set_multi_bit",
}

var scenarioExtNames = map[byte]string{
	0x03: "scenario_menu_ext3",
	0x1d: "scenario_menu_ext1d",
	0x3d: "scenario_menu_ext3d",
}

var op5DNames = map[byte]string{
	0x01: "op_5d_01",
}

var op5FNames = map[byte]string{
	0x01: "op_5f_01",
}

var systemNames = map[byte]string{
	0x02: "load_game",
	0x03: "save_game",
	0x04: "set_title",
	0x05: "make_popup",
	0x20: "game_end",
	0x30: "get_save_title",
	0x31: "check_save_data",
	0x35: "system_unknown1",
	0x36: "system_unknown2",
	0x37: "system_unknown3",
}

var nameNames = map[byte]string{
	0x01: "name_input_box",
	0x02: "name_input_finish",
	0x03: "name_input_start",
	0x04: "name_input_close",
	0x10: "get_name",
	0x11: "set_name",
	0x12: "get_name2",
	0x20: "name_input_dialog",
	0x21: "name_unknown1",
	0x24: "name_input_dialog_multi",
	0x30: "name_unknown2",
	0x31: "name_unknown3",
}

var bufferRegionNames = map[byte]string{
	0x02: "buffer_region_clear_rect",
	0x04: "buffer_region_draw_rect_line",
	0x07: "buffer_region_invert_color",
	0x10: "buffer_region_color_mask",
	0x11: "buffer_region_fadeout_color",
	0x12: "buffer_region_fadeout_color2",
	0x15: "buffer_region_fadeout_color3",
	0x20: "buffer_region_make_mono",
	0x30: "buffer_region_stretch_blit",
	0x32: "buffer_region_stretch_blit_effect",
}

var bufferNames = map[byte]string{
	0x00: "buffer_copy_same_pos",
	0x01: "buffer_copy_new_pos",
	0x02: "buffer_copy_new_pos_mask",
	0x03: "buffer_copy_color",
	0x05: "buffer_swap",
	0x08: "buffer_copy_with_mask",
	0x11: "buffer_copy_whole_screen",
	0x12: "buffer_copy_whole_screen_mask",
	0x20: "buffer_display_strings",
	0x21: "buffer_display_strings_mask",
	0x22: "buffer_display_strings_color",
}

var flashNames = map[byte]string{
	0x01: "flash_fill_color",
	0x10: "flash_screen",
}

var multiPdtNames = map[byte]string{
	0x03: "multi_pdt_slideshow",
	0x04: "multi_pdt_slideshow_loop",
	0x05: "multi_pdt_stop_slideshow_loop",
	0x10: "multi_pdt_scroll",
	0x20: "multi_pdt_scroll2",
	0x30: "multi_pdt_scroll_cancelable",
}

var areaNames = map[byte]string{
	0x02: "area_read_cur_ard",
	0x03: "area_init",
	0x04: "area_get_clicked",
	0x05: "area_get_clicked2",
	0x10: "area_disable",
	0x11: "area_enable",
	0x15: "area_get",
	0x20: "area_assign",
}

var mouseNames = map[byte]string{
	0x01: "mouse_wait_click",
	0x02: "mouse_set_pos",
	0x03: "mouse_flush_click",
	0x20: "cursor_off",
	0x21: "cursor_on",
}

var windowVarNames = map[byte]string{
	0x01: "get_bg_flag_color",
	0x02: "set_bg_flag_color",
	0x03: "get_window_move",
	0x04: "set_window_move",
	0x05: "get_window_clear_box",
	0x06: "set_window_clear_box",
	0x10: "get_window_waku",
	0x11: "set_window_waku",
}

var messageWinNames = map[byte]string{
	0x01: "get_window_msg_pos",
	0x02: "get_window_com_pos",
	0x03: "get_window_sys_pos",
	0x04: "get_window_sub_pos",
	0x05: "get_window_grp_pos",
	0x11: "set_window_msg_pos",
	0x12: "set_window_com_pos",
	0x13: "set_window_sys_pos",
	0x14: "set_window_sub_pos",
	0x15: "set_window_grp_pos",
}

var systemVarNames = map[byte]string{
	0x01: "get_message_size",
	0x02: "set_message_size",
	0x05: "get_msg_moji_size",
	0x06: "set_msg_moji_size",
	0x10: "get_moji_color",
	0x11: "set_moji_color",
	0x12: "get_msg_cancel",
	0x13: "set_msg_cancel",
	0x16: "get_moji_kage",
	0x17: "set_moji_kage",
	0x18: "get_kage_color",
	0x19: "set_kage_color",
	0x1a: "get_sel_cancel",
	0x1b: "set_sel_cancel",
	0x1c: "get_ctrl_key",
	0x1d: "set_ctrl_key",
	0x1e: "get_save_start",
	0x1f: "set_save_start",
	0x20: "get_disable_nvl_text_flag",
	0x21: "set_disable_nvl_text_flag",
	0x22: "get_fade_time",
	0x23: "set_fade_time",
	0x24: "get_cursor_mono",
	0x25: "set_cursor_mono",
	0x26: "get_copy_wind_sw",
	0x27: "set_copy_wind_sw",
	0x28: "get_msg_speed",
	0x29: "set_msg_speed",
	0x2a: "get_msg_speed2",
	0x2b: "set_msg_speed2",
	0x2c: "get_return_key_wait",
	0x2d: "set_return_key_wait",
	0x2e: "get_koe_text_type",
	0x2f: "set_koe_text_type",
	0x30: "get_game_speck_init",
	0x31: "set_cursor_position",
	0x32: "set_disable_key_mouse_flag",
	0x33: "get_game_speck_init2",
	0x34: "set_game_speck_init",
}

var popupNames = map[byte]string{
	0x01: "get_menu_disabled",
	0x02: "set_menu_disabled",
	0x03: "get_item_disabled",
	0x04: "set_item_disabled",
}

var volumeNames = map[byte]string{
	0x01: "get_bgm_volume",
	0x02: "get_wav_volume",
	0x03: "get_koe_volume",
	0x04: "get_se_volume",
	0x11: "set_bgm_volume",
	0x12: "set_wav_volume",
	0x13: "set_koe_volume",
	0x14: "set_se_volume",
	0x21: "mute_bgm",
	0x22: "mute_wav",
	0x23: "mute_koe",
	0x24: "mute_se",
}

var novelNames = map[byte]string{
	0x01: "novel_set_enabled",
	0x02: "novel_unknown1",
	0x03: "novel_unknown2",
	0x04: "novel_unknown3",
	0x05: "novel_unknown4",
	0x10: "novel_value_table",
}

var conditionNames = map[byte]string{
	0x36: "bit_ne",
	0x37: "bit_eq",
	0x38: "ne",
	0x39: "eq",
	0x3a: "flag_ne_const",
	0x3b: "flag_eq_const",
	0x41: "flag_and_const",
	0x42: "flag_and_const2",
	0x43: "flag_xor_const",
	0x44: "flag_gt_const",
	0x45: "flag_lt_const",
	0x46: "flag_ge_const",
	0x47: "flag_le_const",
	0x48: "flag_ne",
	0x49: "flag_eq",
	0x4f: "flag_and",
	0x50: "flag_and2",
	0x51: "flag_xor",
	0x52: "flag_gt",
	0x53: "flag_lt",
	0x54: "flag_ge",
	0x55: "flag_le",
}
